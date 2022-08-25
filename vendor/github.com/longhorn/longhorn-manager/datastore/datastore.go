package datastore

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	batchlisters_v1beta1 "k8s.io/client-go/listers/batch/v1beta1"
	corelisters "k8s.io/client-go/listers/core/v1"
	policylisters "k8s.io/client-go/listers/policy/v1beta1"
	schedulinglisters "k8s.io/client-go/listers/scheduling/v1"
	storagelisters_v1 "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"

	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	lhinformers "github.com/longhorn/longhorn-manager/k8s/pkg/client/informers/externalversions"
	lhlisters "github.com/longhorn/longhorn-manager/k8s/pkg/client/listers/longhorn/v1beta2"
)

var (
	// SkipListerCheck bypass the created longhorn resource validation
	SkipListerCheck = false
)

// DataStore object
type DataStore struct {
	namespace string

	cacheSyncs []cache.InformerSynced

	lhClient                       lhclientset.Interface
	vLister                        lhlisters.VolumeLister
	VolumeInformer                 cache.SharedInformer
	eLister                        lhlisters.EngineLister
	EngineInformer                 cache.SharedInformer
	rLister                        lhlisters.ReplicaLister
	ReplicaInformer                cache.SharedInformer
	iLister                        lhlisters.EngineImageLister
	EngineImageInformer            cache.SharedInformer
	nLister                        lhlisters.NodeLister
	NodeInformer                   cache.SharedInformer
	sLister                        lhlisters.SettingLister
	SettingInformer                cache.SharedInformer
	imLister                       lhlisters.InstanceManagerLister
	InstanceManagerInformer        cache.SharedInformer
	smLister                       lhlisters.ShareManagerLister
	ShareManagerInformer           cache.SharedInformer
	biLister                       lhlisters.BackingImageLister
	BackingImageInformer           cache.SharedInformer
	bimLister                      lhlisters.BackingImageManagerLister
	BackingImageManagerInformer    cache.SharedInformer
	bidsLister                     lhlisters.BackingImageDataSourceLister
	BackingImageDataSourceInformer cache.SharedInformer
	btLister                       lhlisters.BackupTargetLister
	BackupTargetInformer           cache.SharedInformer
	bvLister                       lhlisters.BackupVolumeLister
	BackupVolumeInformer           cache.SharedInformer
	bLister                        lhlisters.BackupLister
	BackupInformer                 cache.SharedInformer
	rjLister                       lhlisters.RecurringJobLister
	RecurringJobInformer           cache.SharedInformer
	oLister                        lhlisters.OrphanLister
	OrphanInformer                 cache.SharedInformer
	snapLister                     lhlisters.SnapshotLister
	SnapshotInformer               cache.SharedInformer

	kubeClient                    clientset.Interface
	pLister                       corelisters.PodLister
	PodInformer                   cache.SharedInformer
	cjLister                      batchlisters_v1beta1.CronJobLister
	CronJobInformer               cache.SharedInformer
	dsLister                      appslisters.DaemonSetLister
	DaemonSetInformer             cache.SharedInformer
	dpLister                      appslisters.DeploymentLister
	DeploymentInformer            cache.SharedInformer
	pvLister                      corelisters.PersistentVolumeLister
	PersistentVolumeInformer      cache.SharedInformer
	pvcLister                     corelisters.PersistentVolumeClaimLister
	PersistentVolumeClaimInformer cache.SharedInformer
	vaLister                      storagelisters_v1.VolumeAttachmentLister
	VolumeAttachmentInformer      cache.SharedInformer
	cfmLister                     corelisters.ConfigMapLister
	ConfigMapInformer             cache.SharedInformer
	secretLister                  corelisters.SecretLister
	SecretInformer                cache.SharedInformer
	knLister                      corelisters.NodeLister
	KubeNodeInformer              cache.SharedInformer
	pcLister                      schedulinglisters.PriorityClassLister
	PriorityClassInformer         cache.SharedInformer
	csiDriverLister               storagelisters_v1.CSIDriverLister
	CSIDriverInformer             cache.SharedInformer
	storageclassLister            storagelisters_v1.StorageClassLister
	StorageClassInformer          cache.SharedInformer
	pdbLister                     policylisters.PodDisruptionBudgetLister
	PodDistrptionBudgetInformer   cache.SharedInformer
	svLister                      corelisters.ServiceLister
	ServiceInformer               cache.SharedInformer
}

// NewDataStore creates new DataStore object
func NewDataStore(
	lhInformerFactory lhinformers.SharedInformerFactory,
	lhClient lhclientset.Interface,
	kubeInformerFactory informers.SharedInformerFactory,
	kubeClient clientset.Interface,
	namespace string) *DataStore {

	cacheSyncs := []cache.InformerSynced{}

	replicaInformer := lhInformerFactory.Longhorn().V1beta2().Replicas()
	cacheSyncs = append(cacheSyncs, replicaInformer.Informer().HasSynced)
	engineInformer := lhInformerFactory.Longhorn().V1beta2().Engines()
	cacheSyncs = append(cacheSyncs, engineInformer.Informer().HasSynced)
	volumeInformer := lhInformerFactory.Longhorn().V1beta2().Volumes()
	cacheSyncs = append(cacheSyncs, volumeInformer.Informer().HasSynced)
	engineImageInformer := lhInformerFactory.Longhorn().V1beta2().EngineImages()
	cacheSyncs = append(cacheSyncs, engineImageInformer.Informer().HasSynced)
	nodeInformer := lhInformerFactory.Longhorn().V1beta2().Nodes()
	cacheSyncs = append(cacheSyncs, nodeInformer.Informer().HasSynced)
	settingInformer := lhInformerFactory.Longhorn().V1beta2().Settings()
	cacheSyncs = append(cacheSyncs, settingInformer.Informer().HasSynced)
	imInformer := lhInformerFactory.Longhorn().V1beta2().InstanceManagers()
	cacheSyncs = append(cacheSyncs, imInformer.Informer().HasSynced)
	smInformer := lhInformerFactory.Longhorn().V1beta2().ShareManagers()
	cacheSyncs = append(cacheSyncs, smInformer.Informer().HasSynced)
	biInformer := lhInformerFactory.Longhorn().V1beta2().BackingImages()
	cacheSyncs = append(cacheSyncs, biInformer.Informer().HasSynced)
	bimInformer := lhInformerFactory.Longhorn().V1beta2().BackingImageManagers()
	cacheSyncs = append(cacheSyncs, bimInformer.Informer().HasSynced)
	bidsInformer := lhInformerFactory.Longhorn().V1beta2().BackingImageDataSources()
	cacheSyncs = append(cacheSyncs, bidsInformer.Informer().HasSynced)
	btInformer := lhInformerFactory.Longhorn().V1beta2().BackupTargets()
	cacheSyncs = append(cacheSyncs, btInformer.Informer().HasSynced)
	bvInformer := lhInformerFactory.Longhorn().V1beta2().BackupVolumes()
	cacheSyncs = append(cacheSyncs, bvInformer.Informer().HasSynced)
	bInformer := lhInformerFactory.Longhorn().V1beta2().Backups()
	cacheSyncs = append(cacheSyncs, bInformer.Informer().HasSynced)
	rjInformer := lhInformerFactory.Longhorn().V1beta2().RecurringJobs()
	cacheSyncs = append(cacheSyncs, rjInformer.Informer().HasSynced)
	oInformer := lhInformerFactory.Longhorn().V1beta2().Orphans()
	cacheSyncs = append(cacheSyncs, oInformer.Informer().HasSynced)
	snapInformer := lhInformerFactory.Longhorn().V1beta2().Snapshots()
	cacheSyncs = append(cacheSyncs, snapInformer.Informer().HasSynced)

	podInformer := kubeInformerFactory.Core().V1().Pods()
	cacheSyncs = append(cacheSyncs, podInformer.Informer().HasSynced)
	kubeNodeInformer := kubeInformerFactory.Core().V1().Nodes()
	cacheSyncs = append(cacheSyncs, kubeNodeInformer.Informer().HasSynced)
	persistentVolumeInformer := kubeInformerFactory.Core().V1().PersistentVolumes()
	cacheSyncs = append(cacheSyncs, persistentVolumeInformer.Informer().HasSynced)
	persistentVolumeClaimInformer := kubeInformerFactory.Core().V1().PersistentVolumeClaims()
	cacheSyncs = append(cacheSyncs, persistentVolumeClaimInformer.Informer().HasSynced)
	volumeAttachmentInformer := kubeInformerFactory.Storage().V1().VolumeAttachments()
	cacheSyncs = append(cacheSyncs, volumeAttachmentInformer.Informer().HasSynced)
	configMapInformer := kubeInformerFactory.Core().V1().ConfigMaps()
	cacheSyncs = append(cacheSyncs, configMapInformer.Informer().HasSynced)
	secretInformer := kubeInformerFactory.Core().V1().Secrets()
	cacheSyncs = append(cacheSyncs, secretInformer.Informer().HasSynced)
	cronJobInformer := kubeInformerFactory.Batch().V1beta1().CronJobs()
	cacheSyncs = append(cacheSyncs, cronJobInformer.Informer().HasSynced)
	daemonSetInformer := kubeInformerFactory.Apps().V1().DaemonSets()
	cacheSyncs = append(cacheSyncs, daemonSetInformer.Informer().HasSynced)
	deploymentInformer := kubeInformerFactory.Apps().V1().Deployments()
	cacheSyncs = append(cacheSyncs, deploymentInformer.Informer().HasSynced)
	priorityClassInformer := kubeInformerFactory.Scheduling().V1().PriorityClasses()
	cacheSyncs = append(cacheSyncs, priorityClassInformer.Informer().HasSynced)
	csiDriverInformer := kubeInformerFactory.Storage().V1().CSIDrivers()
	cacheSyncs = append(cacheSyncs, csiDriverInformer.Informer().HasSynced)
	storageclassInformer := kubeInformerFactory.Storage().V1().StorageClasses()
	cacheSyncs = append(cacheSyncs, storageclassInformer.Informer().HasSynced)
	pdbInformer := kubeInformerFactory.Policy().V1beta1().PodDisruptionBudgets()
	cacheSyncs = append(cacheSyncs, pdbInformer.Informer().HasSynced)
	serviceInformer := kubeInformerFactory.Core().V1().Services()
	cacheSyncs = append(cacheSyncs, serviceInformer.Informer().HasSynced)

	return &DataStore{
		namespace: namespace,

		cacheSyncs: cacheSyncs,

		lhClient:                       lhClient,
		vLister:                        volumeInformer.Lister(),
		VolumeInformer:                 volumeInformer.Informer(),
		eLister:                        engineInformer.Lister(),
		EngineInformer:                 engineInformer.Informer(),
		rLister:                        replicaInformer.Lister(),
		ReplicaInformer:                replicaInformer.Informer(),
		iLister:                        engineImageInformer.Lister(),
		EngineImageInformer:            engineImageInformer.Informer(),
		nLister:                        nodeInformer.Lister(),
		NodeInformer:                   nodeInformer.Informer(),
		sLister:                        settingInformer.Lister(),
		SettingInformer:                settingInformer.Informer(),
		imLister:                       imInformer.Lister(),
		InstanceManagerInformer:        imInformer.Informer(),
		smLister:                       smInformer.Lister(),
		ShareManagerInformer:           smInformer.Informer(),
		biLister:                       biInformer.Lister(),
		BackingImageInformer:           biInformer.Informer(),
		bimLister:                      bimInformer.Lister(),
		BackingImageManagerInformer:    bimInformer.Informer(),
		bidsLister:                     bidsInformer.Lister(),
		BackingImageDataSourceInformer: bidsInformer.Informer(),
		btLister:                       btInformer.Lister(),
		BackupTargetInformer:           btInformer.Informer(),
		bvLister:                       bvInformer.Lister(),
		BackupVolumeInformer:           bvInformer.Informer(),
		bLister:                        bInformer.Lister(),
		BackupInformer:                 bInformer.Informer(),
		rjLister:                       rjInformer.Lister(),
		RecurringJobInformer:           rjInformer.Informer(),
		oLister:                        oInformer.Lister(),
		OrphanInformer:                 oInformer.Informer(),
		snapLister:                     snapInformer.Lister(),
		SnapshotInformer:               snapInformer.Informer(),

		kubeClient:                    kubeClient,
		pLister:                       podInformer.Lister(),
		PodInformer:                   podInformer.Informer(),
		cjLister:                      cronJobInformer.Lister(),
		CronJobInformer:               cronJobInformer.Informer(),
		dsLister:                      daemonSetInformer.Lister(),
		DaemonSetInformer:             daemonSetInformer.Informer(),
		dpLister:                      deploymentInformer.Lister(),
		DeploymentInformer:            deploymentInformer.Informer(),
		pvLister:                      persistentVolumeInformer.Lister(),
		PersistentVolumeInformer:      persistentVolumeInformer.Informer(),
		pvcLister:                     persistentVolumeClaimInformer.Lister(),
		PersistentVolumeClaimInformer: persistentVolumeClaimInformer.Informer(),
		vaLister:                      volumeAttachmentInformer.Lister(),
		VolumeAttachmentInformer:      volumeAttachmentInformer.Informer(),
		cfmLister:                     configMapInformer.Lister(),
		ConfigMapInformer:             configMapInformer.Informer(),
		secretLister:                  secretInformer.Lister(),
		SecretInformer:                secretInformer.Informer(),
		knLister:                      kubeNodeInformer.Lister(),
		KubeNodeInformer:              kubeNodeInformer.Informer(),
		pcLister:                      priorityClassInformer.Lister(),
		PriorityClassInformer:         priorityClassInformer.Informer(),
		csiDriverLister:               csiDriverInformer.Lister(),
		CSIDriverInformer:             csiDriverInformer.Informer(),
		storageclassLister:            storageclassInformer.Lister(),
		StorageClassInformer:          storageclassInformer.Informer(),
		pdbLister:                     pdbInformer.Lister(),
		PodDistrptionBudgetInformer:   pdbInformer.Informer(),
		svLister:                      serviceInformer.Lister(),
		ServiceInformer:               serviceInformer.Informer(),
	}
}

// Sync returns WaitForCacheSync for Longhorn DataStore
func (s *DataStore) Sync(stopCh <-chan struct{}) bool {
	return cache.WaitForNamedCacheSync("longhorn datastore", stopCh, s.cacheSyncs...)
}

// ErrorIsNotFound checks if given error match
// metav1.StatusReasonNotFound
func ErrorIsNotFound(err error) bool {
	return apierrors.IsNotFound(err)
}

// ErrorIsConflict checks if given error match
// metav1.StatusReasonConflict
func ErrorIsConflict(err error) bool {
	return apierrors.IsConflict(err)
}
