package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/upgrade"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	CRDEngineName                 = "engines.longhorn.io"
	CRDReplicaName                = "replicas.longhorn.io"
	CRDVolumeName                 = "volumes.longhorn.io"
	CRDEngineImageName            = "engineimages.longhorn.io"
	CRDNodeName                   = "nodes.longhorn.io"
	CRDInstanceManagerName        = "instancemanagers.longhorn.io"
	CRDShareManagerName           = "sharemanagers.longhorn.io"
	CRDBackingImageName           = "backingimages.longhorn.io"
	CRDBackingImageManagerName    = "backingimagemanagers.longhorn.io"
	CRDBackingImageDataSourceName = "backingimagedatasources.longhorn.io"
	CRDBackupTargetName           = "backuptargets.longhorn.io"
	CRDBackupVolumeName           = "backupvolumes.longhorn.io"
	CRDBackupName                 = "backups.longhorn.io"
	CRDRecurringJobName           = "recurringjobs.longhorn.io"
	CRDOrphanName                 = "orphans.longhorn.io"
	CRDSnapshotName               = "snapshots.longhorn.io"

	EnvLonghornNamespace = "LONGHORN_NAMESPACE"
)

var (
	gracePeriod = 90 * time.Second
)

type UninstallController struct {
	*baseController
	namespace string
	force     bool
	ds        *datastore.DataStore
	stopCh    chan struct{}

	kubeClient clientset.Interface

	cacheSyncs []cache.InformerSynced
}

func NewUninstallController(
	logger logrus.FieldLogger,
	namespace string,
	force bool,
	ds *datastore.DataStore,
	stopCh chan struct{},
	kubeClient clientset.Interface,
	extensionsClient apiextension.Interface,
) *UninstallController {
	c := &UninstallController{
		baseController: newBaseControllerWithQueue("longhorn-uninstall", logger,
			workqueue.NewNamedRateLimitingQueue(workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemExponentialFailureRateLimiter(100*time.Millisecond, 2*time.Second),
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(100), 1000)},
			), "longhorn-uninstall"),
		),
		namespace: namespace,
		force:     force,
		ds:        ds,
		stopCh:    stopCh,

		kubeClient: kubeClient,
	}

	ds.CSIDriverInformer.AddEventHandler(c.controlleeHandler())
	ds.DaemonSetInformer.AddEventHandler(c.namespacedControlleeHandler())
	ds.DeploymentInformer.AddEventHandler(c.namespacedControlleeHandler())
	cacheSyncs := []cache.InformerSynced{
		ds.CSIDriverInformer.HasSynced,
		ds.DaemonSetInformer.HasSynced,
		ds.DeploymentInformer.HasSynced,
	}

	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDEngineName, metav1.GetOptions{}); err == nil {
		ds.EngineInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.EngineInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDReplicaName, metav1.GetOptions{}); err == nil {
		ds.ReplicaInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.ReplicaInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDVolumeName, metav1.GetOptions{}); err == nil {
		ds.VolumeInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.VolumeInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDEngineImageName, metav1.GetOptions{}); err == nil {
		ds.EngineImageInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.EngineImageInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDNodeName, metav1.GetOptions{}); err == nil {
		ds.NodeInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.NodeInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDInstanceManagerName, metav1.GetOptions{}); err == nil {
		ds.InstanceManagerInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.InstanceManagerInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDShareManagerName, metav1.GetOptions{}); err == nil {
		ds.ShareManagerInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.ShareManagerInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDBackingImageName, metav1.GetOptions{}); err == nil {
		ds.BackingImageInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.BackingImageInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDBackingImageManagerName, metav1.GetOptions{}); err == nil {
		ds.BackingImageManagerInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.BackingImageManagerInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDBackingImageDataSourceName, metav1.GetOptions{}); err == nil {
		ds.BackingImageDataSourceInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.BackingImageDataSourceInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDBackupTargetName, metav1.GetOptions{}); err == nil {
		ds.BackupTargetInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.BackupTargetInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDBackupVolumeName, metav1.GetOptions{}); err == nil {
		ds.BackupVolumeInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.BackupVolumeInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDBackupName, metav1.GetOptions{}); err == nil {
		ds.BackupInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.BackupInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDRecurringJobName, metav1.GetOptions{}); err == nil {
		ds.RecurringJobInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.RecurringJobInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDOrphanName, metav1.GetOptions{}); err == nil {
		ds.OrphanInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.OrphanInformer.HasSynced)
	}
	if _, err := extensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), CRDSnapshotName, metav1.GetOptions{}); err == nil {
		ds.SnapshotInformer.AddEventHandler(c.controlleeHandler())
		cacheSyncs = append(cacheSyncs, ds.SnapshotInformer.HasSynced)
	}

	c.cacheSyncs = cacheSyncs

	return c
}

func (c *UninstallController) controlleeHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.enqueueControlleeChange() },
		UpdateFunc: func(old, cur interface{}) { c.enqueueControlleeChange() },
		DeleteFunc: func(obj interface{}) { c.enqueueControlleeChange() },
	}
}

func (c *UninstallController) namespacedControlleeHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.enqueueNamespacedControlleeChange(obj) },
		UpdateFunc: func(old, cur interface{}) { c.enqueueNamespacedControlleeChange(cur) },
		DeleteFunc: func(obj interface{}) { c.enqueueNamespacedControlleeChange(obj) },
	}
}

func (c *UninstallController) enqueueControlleeChange() {
	c.queue.Add("uninstall")
}

func (c *UninstallController) enqueueNamespacedControlleeChange(obj interface{}) {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get meta for object %#v: %v", obj, err))
		return
	}

	if metadata.GetNamespace() == os.Getenv(EnvLonghornNamespace) {
		c.enqueueControlleeChange()
	}
}

func (c *UninstallController) Run() error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	if !cache.WaitForNamedCacheSync("longhorn uninstall", c.stopCh, c.cacheSyncs...) {
		return fmt.Errorf("failed to sync informers")
	}

	if err := c.checkPreconditions(); err != nil {
		close(c.stopCh)
		return err
	}

	startTime := time.Now()
	c.logger.Info("Uninstalling...")
	defer func() {
		log := c.logger.WithField("runtime", time.Since(startTime))
		log.Info("Uninstallation completed")
	}()
	go wait.Until(c.worker, time.Second, c.stopCh)

	<-c.stopCh
	return nil
}

func (c *UninstallController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *UninstallController) processNextWorkItem() bool {
	key, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.uninstall()
	c.handleErr(err, key)

	return true
}

func (c *UninstallController) handleErr(err error, key interface{}) {
	if err == nil {
		c.queue.Forget(key)
		return
	}

	c.logger.WithError(err).Warn("worker error")
	c.queue.AddRateLimited(key)
}

func getLoggerForUninstallCSIDriver(logger logrus.FieldLogger, name string) *logrus.Entry {
	return logger.WithField("CSIDriver", name)
}

func getLoggerForUninstallDaemonSet(logger logrus.FieldLogger, name string) *logrus.Entry {
	return logger.WithField("daemonSet", name)
}

func getLoggerForUninstallDeployment(logger logrus.FieldLogger, name string) *logrus.Entry {
	return logger.WithField("deployment", name)
}

func getLoggerForUninstallService(logger logrus.FieldLogger, name string) *logrus.Entry {
	return logger.WithField("service", name)
}

func (c *UninstallController) uninstall() error {
	if ready, err := c.managerReady(); err != nil {
		return err
	} else if ready {
		if waitForUpdate, err := c.deleteCRDs(); err != nil || waitForUpdate {
			return err
		}
	}

	// A race condition exists where manager may attempt to recreate certain
	// CRDs after their deletion. We must delete manager and do a final check
	// for CRDs, just in case.
	if waitForUpdate, err := c.deleteManager(); err != nil || waitForUpdate {
		return err
	}

	if waitForUpdate, err := c.deleteDriver(); err != nil || waitForUpdate {
		return err
	}

	// We set gracePeriod=0s because there is no possibility of graceful
	// cleanup without a running manager.
	gracePeriod = 0 * time.Second
	if waitForUpdate, err := c.deleteCRDs(); err != nil || waitForUpdate {
		return err
	}

	if waitForUpdate, err := c.deleteWebhookDeployment(types.LonghornAdmissionWebhookDeploymentName); err != nil || waitForUpdate {
		return err
	}

	if waitForUpdate, err := c.deleteWebhookDeployment(types.LonghornConversionWebhookDeploymentName); err != nil || waitForUpdate {
		return err
	}

	if err := c.deleteWebhookConfiguration(); err != nil {
		return err
	}

	if err := c.deleteStorageClass(); err != nil {
		return err
	}

	if err := c.deleteLease(); err != nil {
		return err
	}

	// Success
	close(c.stopCh)
	return nil
}

func (c *UninstallController) checkPreconditions() error {
	if ready, err := c.managerReady(); err != nil {
		return err
	} else if !ready {
		if c.force {
			c.logger.Warn("Manager not ready, this may leave data behind")
			gracePeriod = 0 * time.Second
		} else {
			return fmt.Errorf("manager not ready, set --force to continue")
		}
	}

	if vols, err := c.ds.ListVolumesRO(); err != nil {
		return err
	} else if len(vols) > 0 {
		volumesInUse := false
		for _, vol := range vols {
			if vol.Status.State == longhorn.VolumeStateAttaching ||
				vol.Status.State == longhorn.VolumeStateAttached {
				log := getLoggerForVolume(c.logger, vol)
				log.Warn("Volume in use")
				volumesInUse = true
			}
		}
		if volumesInUse && !c.force {
			return fmt.Errorf("volume(s) in use, set --force to continue")
		}
	}

	return nil
}

func (c *UninstallController) deleteStorageClass() error {
	err := c.ds.DeleteStorageClass(types.DefaultStorageClassName)
	if err != nil && !datastore.ErrorIsNotFound(err) {
		return err
	}
	return nil
}

func (c *UninstallController) deleteLease() error {
	err := c.ds.DeleteLease(upgrade.LeaseLockName)
	if err != nil && !datastore.ErrorIsNotFound(err) {
		return err
	}
	return nil
}

func (c *UninstallController) deleteCRDs() (bool, error) {
	if volumes, err := c.ds.ListVolumes(); err != nil {
		return true, err
	} else if len(volumes) > 0 {
		c.logger.Infof("Found %d volumes remaining", len(volumes))
		return true, c.deleteVolumes(volumes)
	}

	if snapshots, err := c.ds.ListSnapshots(); err != nil {
		return true, err
	} else if len(snapshots) > 0 {
		// We deleted all volume CRs before deleting snapshot CRs in the above steps.
		// Since at this step the volume is already gone, we can delete all snapshot CRs in the system
		c.logger.Infof("Found %d snapshots remaining", len(snapshots))
		return true, c.deleteSnapshots(snapshots)
	}

	if engines, err := c.ds.ListEngines(); err != nil {
		return true, err
	} else if len(engines) > 0 {
		c.logger.Infof("Found %d engines remaining", len(engines))
		return true, c.deleteEngines(engines)
	}

	if replicas, err := c.ds.ListReplicas(); err != nil {
		return true, err
	} else if len(replicas) > 0 {
		c.logger.Infof("Found %d replicas remaining", len(replicas))
		return true, c.deleteReplicas(replicas)
	}

	// Unset backup target to prevent the remote backup target
	// backup volume config, and backup config and it's data
	// being deleted during uninstall process.
	// TODO:
	//   Remove the setting CRs and add OwnerReferences on BackupTarget CR.
	//   After that when deleting setting CRs, the default BackupTarget CR
	//   be cascading deleted automatically.
	targetSetting, err := c.ds.GetSetting(types.SettingNameBackupTarget)
	if err != nil {
		return true, err
	}
	if targetSetting.Value != "" {
		targetSetting.Value = ""
		if _, err := c.ds.UpdateSetting(targetSetting); err != nil {
			return true, err
		}
	}

	// Waits the BackupVolume CRs be clean up by backup_target_controller
	if backupVolumes, err := c.ds.ListBackupVolumes(); err != nil {
		return true, err
	} else if len(backupVolumes) > 0 {
		c.logger.Infof("Found %d backupvolumes remaining", len(backupVolumes))
		return true, nil
	}

	// Waits the Backup CRs be clean up by backup_volume_controller
	if backups, err := c.ds.ListBackups(); err != nil {
		return true, err
	} else if len(backups) > 0 {
		c.logger.Infof("Found %d backups remaining", len(backups))
		return true, nil
	}

	// Delete the BackupTarget CRs
	if backupTargets, err := c.ds.ListBackupTargets(); err != nil {
		return true, err
	} else if len(backupTargets) > 0 {
		c.logger.Infof("Found %d backuptargets remaining", len(backupTargets))
		return true, c.deleteBackupTargets(backupTargets)
	}

	if engineImages, err := c.ds.ListEngineImages(); err != nil {
		return true, err
	} else if len(engineImages) > 0 {
		c.logger.Infof("Found %d engineimages remaining", len(engineImages))
		return true, c.deleteEngineImages(engineImages)
	}

	if backingImages, err := c.ds.ListBackingImages(); err != nil {
		return true, err
	} else if len(backingImages) > 0 {
		c.logger.Infof("Found %d backingimages remaining", len(backingImages))
		return true, c.deleteBackingImages(backingImages)
	}

	if shareManagers, err := c.ds.ListShareManagers(); err != nil {
		return true, err
	} else if len(shareManagers) > 0 {
		c.logger.Infof("Found %d share managers remaining", len(shareManagers))
		return true, c.deleteShareManagers(shareManagers)
	}

	if backingImageManagers, err := c.ds.ListBackingImageManagers(); err != nil {
		return true, err
	} else if len(backingImageManagers) > 0 {
		c.logger.Infof("Found %d backingimagemanagers remaining", len(backingImageManagers))
		return true, c.deleteBackingImageManagers(backingImageManagers)
	}

	if backingImageDataSources, err := c.ds.ListBackingImageDataSources(); err != nil {
		return true, err
	} else if len(backingImageDataSources) > 0 {
		c.logger.Infof("Found %d backingImageDataSources remaining", len(backingImageDataSources))
		return true, c.deleteBackingImageDataSource(backingImageDataSources)
	}

	if recurringJobs, err := c.ds.ListRecurringJobs(); err != nil {
		return true, err
	} else if len(recurringJobs) > 0 {
		c.logger.Infof("Found %d recurring jobs remaining", len(recurringJobs))
		return true, c.deleteRecurringJobs(recurringJobs)
	}

	if nodes, err := c.ds.ListNodes(); err != nil {
		return true, err
	} else if len(nodes) > 0 {
		c.logger.Infof("Found %d nodes remaining", len(nodes))
		return true, c.deleteNodes(nodes)
	}

	if instanceManagers, err := c.ds.ListInstanceManagers(); err != nil {
		return true, err
	} else if len(instanceManagers) > 0 {
		c.logger.Infof("Found %d instance managers remaining", len(instanceManagers))
		return true, c.deleteInstanceManagers(instanceManagers)
	}

	if orphans, err := c.ds.ListOrphans(); err != nil {
		return true, err
	} else if len(orphans) > 0 {
		c.logger.Infof("Found %d orphans remaining", len(orphans))
		return true, c.deleteOrphans(orphans)
	}

	return false, nil
}

func (c *UninstallController) deleteVolumes(vols map[string]*longhorn.Volume) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete volumes")
	}()
	for _, vol := range vols {
		log := getLoggerForVolume(c.logger, vol)

		timeout := metav1.NewTime(time.Now().Add(-gracePeriod))
		if vol.DeletionTimestamp == nil {
			if err = c.ds.DeleteVolume(vol.Name); err != nil {
				err = errors.Wrapf(err, "Failed to mark for deletion")
				return
			}
			log.Info("Marked for deletion")
		} else if vol.DeletionTimestamp.Before(&timeout) {
			if err = c.ds.RemoveFinalizerForVolume(vol); err != nil {
				err = errors.Wrapf(err, "Failed to remove finalizer")
				return
			}
			log.Info("Removed finalizer")
		}
	}
	return
}

func (c *UninstallController) deleteSnapshots(snapshots map[string]*longhorn.Snapshot) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete snapshots")
	}()
	for _, snap := range snapshots {
		log := getLoggerForSnapshot(c.logger, snap)

		timeout := metav1.NewTime(time.Now().Add(-gracePeriod))
		if snap.DeletionTimestamp == nil {
			if err = c.ds.DeleteSnapshot(snap.Name); err != nil {
				err = errors.Wrapf(err, "Failed to mark for deletion")
				return
			}
			log.Info("Marked for deletion")
		} else if snap.DeletionTimestamp.Before(&timeout) {
			if err = c.ds.RemoveFinalizerForSnapshot(snap); err != nil {
				err = errors.Wrapf(err, "Failed to remove finalizer")
				return
			}
			log.Info("Removed finalizer")
		}
	}
	return
}

func (c *UninstallController) deleteEngines(engines map[string]*longhorn.Engine) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete engines")
	}()
	for _, engine := range engines {
		log := getLoggerForEngine(c.logger, engine)

		timeout := metav1.NewTime(time.Now().Add(-gracePeriod))
		if engine.DeletionTimestamp == nil {
			if err = c.ds.DeleteEngine(engine.Name); err != nil {
				err = errors.Wrapf(err, "Failed to mark for deletion")
				return
			}
			log.Info("Marked for deletion")
		} else if engine.DeletionTimestamp.Before(&timeout) {
			if err = c.ds.RemoveFinalizerForEngine(engine); err != nil {
				err = errors.Wrapf(err, "Failed to remove finalizer")
				return
			}
			log.Info("Removed finalizer")
		}
	}
	return
}

func (c *UninstallController) deleteReplicas(replicas map[string]*longhorn.Replica) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete replicas")
	}()
	for _, replica := range replicas {
		log := getLoggerForReplica(c.logger, replica)

		timeout := metav1.NewTime(time.Now().Add(-gracePeriod))
		if replica.DeletionTimestamp == nil {
			if err = c.ds.DeleteReplica(replica.Name); err != nil {
				err = errors.Wrapf(err, "Failed to mark for deletion")
				return
			}
			log.Info("Marked for deletion")
		} else if replica.DeletionTimestamp.Before(&timeout) {
			if err = c.ds.RemoveFinalizerForReplica(replica); err != nil {
				err = errors.Wrapf(err, "Failed to remove finalizer")
				return
			}
			log.Info("Removed finalizer")
		}
	}
	return
}

func (c *UninstallController) deleteBackupTargets(backupTargets map[string]*longhorn.BackupTarget) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete backup targets")
	}()
	for _, bt := range backupTargets {
		log := getLoggerForBackupTarget(c.logger, bt)
		if bt.DeletionTimestamp == nil {
			if err = c.ds.DeleteBackupTarget(bt.Name); err != nil {
				return errors.Wrapf(err, "Failed to mark for deletion")
			}
			log.Info("Marked for deletion")
		}
	}
	return
}

func (c *UninstallController) deleteEngineImages(engineImages map[string]*longhorn.EngineImage) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete engine images")
	}()
	for _, ei := range engineImages {
		log := getLoggerForEngineImage(c.logger, ei)

		timeout := metav1.NewTime(time.Now().Add(-gracePeriod))
		if ei.DeletionTimestamp == nil {
			if err = c.ds.DeleteEngineImage(ei.Name); err != nil {
				err = errors.Wrapf(err, "Failed to mark for deletion")
				return
			}
			log.Info("Marked for deletion")
		} else if ei.DeletionTimestamp.Before(&timeout) {
			dsName := types.GetDaemonSetNameFromEngineImageName(ei.Name)
			if err = c.ds.DeleteDaemonSet(dsName); err != nil {
				if !apierrors.IsNotFound(err) {
					err = errors.Wrapf(err, "Failed to remove daemon set")
					return
				}
				log.Info("Removed daemon set")
				err = nil
			}
			if err = c.ds.RemoveFinalizerForEngineImage(ei); err != nil {
				err = errors.Wrapf(err, "Failed to remove finalizer")
				return
			}
			log.Info("Removed finalizer")
		}
	}
	return
}

func (c *UninstallController) deleteNodes(nodes map[string]*longhorn.Node) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete nodes")
	}()
	for _, node := range nodes {
		log := getLoggerForNode(c.logger, node)

		if node.DeletionTimestamp == nil {
			if err = c.ds.DeleteNode(node.Name); err != nil {
				err = errors.Wrapf(err, "Failed to mark for deletion")
				return
			}
			log.Info("Marked for deletion")
		} else {
			if err = c.ds.RemoveFinalizerForNode(node); err != nil {
				err = errors.Wrapf(err, "Failed to remove finalizer")
				return
			}
			log.Info("Removed finalizer")
		}
	}
	return
}

func (c *UninstallController) deleteInstanceManagers(instanceManagers map[string]*longhorn.InstanceManager) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete instance managers")
	}()
	for _, im := range instanceManagers {
		log := getLoggerForInstanceManager(c.logger, im)
		if im.DeletionTimestamp == nil {
			if err = c.ds.DeleteInstanceManager(im.Name); err != nil {
				err = errors.Wrapf(err, "Failed to mark for deletion")
				return
			}
			log.Info("Marked for deletion")
		}
	}
	return
}

func (c *UninstallController) deleteShareManagers(shareManagers map[string]*longhorn.ShareManager) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete share managers")
	}()
	for _, sm := range shareManagers {
		log := getLoggerForShareManager(c.logger, sm)

		timeout := metav1.NewTime(time.Now().Add(-gracePeriod))
		if sm.DeletionTimestamp == nil {
			if err = c.ds.DeleteShareManager(sm.Name); err != nil {
				err = errors.Wrapf(err, "Failed to mark for deletion")
				return
			}
			log.Info("Marked for deletion")
		} else if sm.DeletionTimestamp.Before(&timeout) {
			podName := types.GetShareManagerPodNameFromShareManagerName(sm.Name)
			if err = c.ds.DeletePod(podName); err != nil {
				return err
			}
			log.Infof("Removing share manager related pod %v", podName)

			if err = c.ds.RemoveFinalizerForShareManager(sm); err != nil {
				err = errors.Wrapf(err, "Failed to remove finalizer")
				return
			}
			log.Info("Removed finalizer")
		}
	}
	return
}

func (c *UninstallController) deleteBackingImages(backingImages map[string]*longhorn.BackingImage) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete backing images")
	}()
	for _, bi := range backingImages {
		log := getLoggerForBackingImage(c.logger, bi)

		timeout := metav1.NewTime(time.Now().Add(-gracePeriod))
		if bi.DeletionTimestamp == nil {
			if err = c.ds.DeleteBackingImage(bi.Name); err != nil {
				return errors.Wrapf(err, "Failed to mark for deletion")
			}
			log.Info("Marked for deletion")
		} else if bi.DeletionTimestamp.Before(&timeout) {
			if err = c.ds.RemoveFinalizerForBackingImage(bi); err != nil {
				return errors.Wrapf(err, "Failed to remove finalizer")
			}
			log.Info("Removed finalizer")
		}
	}
	return nil
}

func (c *UninstallController) deleteBackingImageManagers(backingImageManagers map[string]*longhorn.BackingImageManager) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete backing image managers")
	}()
	for _, bim := range backingImageManagers {
		log := getLoggerForBackingImageManager(c.logger, bim)

		timeout := metav1.NewTime(time.Now().Add(-gracePeriod))
		if bim.DeletionTimestamp == nil {
			if err = c.ds.DeleteBackingImageManager(bim.Name); err != nil {
				return errors.Wrapf(err, "Failed to mark for deletion")
			}
			log.Info("Marked for deletion")
		} else if bim.DeletionTimestamp.Before(&timeout) {
			if err := c.ds.DeletePod(bim.Name); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			if err = c.ds.RemoveFinalizerForBackingImageManager(bim); err != nil {
				return errors.Wrapf(err, "Failed to remove finalizer")
			}
			log.Info("Removed finalizer")
		}
	}
	return nil
}

func (c *UninstallController) deleteBackingImageDataSource(backingImageDataSources map[string]*longhorn.BackingImageDataSource) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete backing image data sources")
	}()
	for _, bids := range backingImageDataSources {
		log := getLoggerForBackingImageDataSource(c.logger, bids)

		timeout := metav1.NewTime(time.Now().Add(-gracePeriod))
		if bids.DeletionTimestamp == nil {
			if err = c.ds.DeleteBackingImageDataSource(bids.Name); err != nil {
				return errors.Wrapf(err, "Failed to mark for deletion")
			}
			log.Info("Marked for deletion")
		} else if bids.DeletionTimestamp.Before(&timeout) {
			if err := c.ds.DeletePod(bids.Name); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			if err = c.ds.RemoveFinalizerForBackingImageDataSource(bids); err != nil {
				return errors.Wrapf(err, "Failed to remove finalizer")
			}
			log.Info("Removed finalizer")
		}
	}
	return nil
}

func (c *UninstallController) deleteRecurringJobs(recurringJobs map[string]*longhorn.RecurringJob) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete recurring jobs")
	}()
	for _, job := range recurringJobs {
		log := getLoggerForRecurringJob(c.logger, job)
		if job.DeletionTimestamp == nil {
			if err = c.ds.DeleteRecurringJob(job.Name); err != nil {
				return errors.Wrapf(err, "Failed to mark for deletion")
			}
			log.Info("Marked for deletion")
		}
	}
	return nil
}

func (c *UninstallController) deleteOrphans(orphans map[string]*longhorn.Orphan) (err error) {
	defer func() {
		err = errors.Wrapf(err, "Failed to delete orphans")
	}()
	for _, orphan := range orphans {
		log := getLoggerForOrphan(c.logger, orphan)
		if orphan.DeletionTimestamp == nil {
			if err = c.ds.DeleteOrphan(orphan.Name); err != nil {
				return errors.Wrapf(err, "Failed to mark for deletion")
			}
			log.Info("Marked for deletion")
		}
	}
	return nil
}

func (c *UninstallController) deleteManager() (bool, error) {
	log := getLoggerForUninstallDaemonSet(c.logger, types.LonghornManagerDaemonSetName)

	if ds, err := c.ds.GetDaemonSet(types.LonghornManagerDaemonSetName); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return true, err
	} else if ds.DeletionTimestamp == nil {
		if err := c.ds.DeleteDaemonSet(types.LonghornManagerDaemonSetName); err != nil {
			log.Warn("failed to mark for deletion")
			return true, err
		}
		log.Info("Marked for deletion")
		return true, nil
	}
	log.Info("Already marked for deletion")
	return true, nil
}

func (c *UninstallController) deleteWebhookDeployment(deployment string) (bool, error) {
	log := getLoggerForUninstallDeployment(c.logger, deployment)

	if ds, err := c.ds.GetDeployment(deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return true, err
	} else if ds.DeletionTimestamp == nil {
		if err := c.ds.DeleteDeployment(deployment); err != nil {
			log.Warn("failed to mark for deletion")
			return true, err
		}
		log.Info("Marked for deletion")
		return true, nil
	}
	log.Info("Already marked for deletion")
	return true, nil
}

func (c *UninstallController) deleteWebhookConfiguration() error {
	validatingCfg, err := c.kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(context.TODO(), types.ValidatingWebhookName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if validatingCfg != nil {
		if err := c.kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(context.TODO(), types.ValidatingWebhookName, metav1.DeleteOptions{}); err != nil {
			return err
		}
		c.logger.Infof("Successfully clean up the validating webhook configuration %s", types.ValidatingWebhookName)
	}

	mutatingCfg, err := c.kubeClient.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.TODO(), types.MutatingWebhookName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if mutatingCfg != nil {
		if err := c.kubeClient.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(context.TODO(), types.MutatingWebhookName, metav1.DeleteOptions{}); err != nil {
			return err
		}
		c.logger.Infof("Successfully clean up the mutating webhook configuration %s", types.MutatingWebhookName)
	}
	return nil
}

func (c *UninstallController) managerReady() (bool, error) {
	log := getLoggerForUninstallDaemonSet(c.logger, types.LonghornManagerDaemonSetName)

	if ds, err := c.ds.GetDaemonSet(types.LonghornManagerDaemonSetName); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	} else if ds.DeletionTimestamp != nil {
		log.Warn("Marked for deletion")
		return false, nil
	} else if ds.Status.NumberReady < ds.Status.DesiredNumberScheduled-1 {
		// During upgrade, there may be at most one pod missing, so we
		// will allow that to support uninstallation during upgrade
		log.Warnf("Not enough ready pods (%d/%d)",
			ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
		return false, nil
	}
	return true, nil
}

func (c *UninstallController) deleteDriver() (bool, error) {
	deploymentsToClean := []string{
		types.DriverDeployerName,
		types.CSIAttacherName,
		types.CSIProvisionerName,
		types.CSIResizerName,
		types.CSISnapshotterName,
	}
	wait := false
	for _, name := range deploymentsToClean {
		log := getLoggerForUninstallDeployment(c.logger, name)

		if driver, err := c.ds.GetDeployment(name); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			log.WithError(err).Warn("Failed to get for deletion")
			wait = true
			continue
		} else if driver.DeletionTimestamp == nil {
			if err := c.ds.DeleteDeployment(name); err != nil {
				log.Warn("Failed to mark for deletion")
				wait = true
				continue
			}
			log.Info("Marked for deletion")
			wait = true
			continue
		}
		log.Info("Already marked for deletion")
		wait = true
	}

	servicesToClean := []string{
		types.CSIAttacherName,
		types.CSIProvisionerName,
		types.CSIResizerName,
		types.CSISnapshotterName,
	}
	for _, name := range servicesToClean {
		log := getLoggerForUninstallService(c.logger, name)

		if service, err := c.ds.GetService(c.namespace, name); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			log.WithError(err).Warn("Failed to get for deletion")
			wait = true
			continue
		} else if service.DeletionTimestamp == nil {
			if err := c.ds.DeleteService(c.namespace, name); err != nil {
				log.Warn("Failed to mark for deletion")
				wait = true
				continue
			}
			log.Info("Marked for deletion")
			wait = true
			continue
		}
		log.Info("Already marked for deletion")
		wait = true
	}

	daemonSetsToClean := []string{
		types.CSIPluginName,
	}
	for _, name := range daemonSetsToClean {
		log := getLoggerForUninstallDaemonSet(c.logger, name)

		if driver, err := c.ds.GetDaemonSet(name); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			log.WithError(err).Warn("Failed to get for deletion")
			wait = true
			continue
		} else if driver.DeletionTimestamp == nil {
			if err := c.ds.DeleteDaemonSet(name); err != nil {
				log.WithError(err).Warn("Failed to mark for deletion")
				wait = true
				continue
			}
			log.Info("Marked for deletion")
			wait = true
			continue
		}
		log.Info("Already marked for deletion")
		wait = true
	}

	if err := c.ds.DeleteCSIDriver(types.LonghornDriverName); err != nil {
		if !apierrors.IsNotFound(err) {
			log := getLoggerForUninstallCSIDriver(c.logger, types.LonghornDriverName)
			log.WithError(err).Warn("Failed to delete")
			wait = true
		}
	}

	return wait, nil
}
