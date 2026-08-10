package imagevolumesizecheck

import (
	"context"
	"fmt"
	"time"

	"github.com/rancher/wrangler/v3/pkg/generated/controllers/core"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/generated/controllers/storage"
	ctlstoragev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/storage/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	ctlharvester "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	ctlkv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/util"
	indexeresutil "github.com/harvester/harvester/pkg/util/indexeres"
	webhookutil "github.com/harvester/harvester/pkg/webhook/util"
)

const (
	imageVolumeSizeCheckCompleted = "ImageVolumeSizeCheckCompleted"
)

// Options configures a Checker.
type Options struct {
	KubeConfigPath string
	KubeContext    string
	Upgrade        string
}

// Checker scans all PVCs cluster-wide for volumes created from a
// VirtualMachineImage whose size is smaller than the image's virtual size,
// and reports them. It is read-only: PVCs can never be shrunk, and
// expanding an in-use volume is a decision best left to the cluster
// operator, so the checker never modifies any resource, it only detects
// and reports violations.
type Checker struct {
	kubeConfig  string
	kubeContext string
	upgradeName string

	harvFactory    *ctlharvester.Factory
	coreFactory    *core.Factory
	virtFactory    *kubevirt.Factory
	storageFactory *storage.Factory

	imageCache    ctlharvesterv1.VirtualMachineImageCache
	upgradeCache  ctlharvesterv1.UpgradeCache
	settingCache  ctlharvesterv1.SettingCache
	pvcCache      ctlcorev1.PersistentVolumeClaimCache
	vmCache       ctlkv1.VirtualMachineCache
	kubevirtCache ctlkv1.KubeVirtCache
	scCache       ctlstoragev1.StorageClassCache

	recorder record.EventRecorder
}

// Assessment summarizes the outcome of a single Run.
type Assessment struct {
	Scanned    int
	Violations []string
}

func NewChecker(options Options) *Checker {
	return &Checker{
		kubeConfig:  options.KubeConfigPath,
		kubeContext: options.KubeContext,
		upgradeName: options.Upgrade,
	}
}

func (c *Checker) Init() error {
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{
			ExplicitPath: c.kubeConfig,
		},
		&clientcmd.ConfigOverrides{
			CurrentContext: c.kubeContext,
		},
	)
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to build REST config: %w", err)
	}

	k8sClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	harvFactory, err := ctlharvester.NewFactoryFromConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create harvester factory: %w", err)
	}
	c.harvFactory = harvFactory
	c.imageCache = harvFactory.Harvesterhci().V1beta1().VirtualMachineImage().Cache()
	c.upgradeCache = harvFactory.Harvesterhci().V1beta1().Upgrade().Cache()
	c.settingCache = harvFactory.Harvesterhci().V1beta1().Setting().Cache()

	coreFactory, err := core.NewFactoryFromConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create core factory: %w", err)
	}
	c.coreFactory = coreFactory
	c.pvcCache = coreFactory.Core().V1().PersistentVolumeClaim().Cache()

	virtFactory, err := kubevirt.NewFactoryFromConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create kubevirt factory: %w", err)
	}
	c.virtFactory = virtFactory
	c.vmCache = virtFactory.Kubevirt().V1().VirtualMachine().Cache()
	c.kubevirtCache = virtFactory.Kubevirt().V1().KubeVirt().Cache()
	// webhookutil.CheckExpand looks up VMs by PVC via this index; it must be
	// registered before the informer starts (see pkg/webhook/indexeres).
	c.vmCache.AddIndexer(indexeresutil.VMByPVCIndex, indexeresutil.VMByPVC)

	storageFactory, err := storage.NewFactoryFromConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create storage factory: %w", err)
	}
	c.storageFactory = storageFactory
	c.scCache = storageFactory.Storage().V1().StorageClass().Cache()

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: k8sClient.CoreV1().Events(util.HarvesterSystemNamespaceName)})
	c.recorder = broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "image-volume-size-check"})

	return nil
}

func (c *Checker) Run(ctx context.Context) (*Assessment, error) {
	defer func() {
		// Wait for events to be flushed
		time.Sleep(10 * time.Second)
	}()

	if err := c.harvFactory.Sync(ctx); err != nil {
		return nil, fmt.Errorf("failed to sync Harvester factory: %w", err)
	}
	if err := c.coreFactory.Sync(ctx); err != nil {
		return nil, fmt.Errorf("failed to sync core factory: %w", err)
	}
	if err := c.virtFactory.Sync(ctx); err != nil {
		return nil, fmt.Errorf("failed to sync KubeVirt factory: %w", err)
	}
	if err := c.storageFactory.Sync(ctx); err != nil {
		return nil, fmt.Errorf("failed to sync storage factory: %w", err)
	}

	pvcs, err := c.pvcCache.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("failed to list PVCs: %w", err)
	}

	assessment := &Assessment{Scanned: len(pvcs)}
	for _, pvc := range pvcs {
		violation, minSize, err := c.checkPVCMinSize(pvc)
		if err != nil {
			logrus.Warnf("failed to check PVC %q: %v", util.GetNamespacedName(pvc), err)
			continue
		}
		if !violation {
			continue
		}

		detail := fmt.Sprintf("%q: current size is smaller than the required minimum of %s", util.GetNamespacedName(pvc), minSize.String())
		if err := webhookutil.CheckExpand(pvc, c.vmCache, c.kubevirtCache, c.scCache, c.settingCache); err != nil {
			detail += fmt.Sprintf("; not expandable right now: %v", err)
		} else {
			detail += "; can be expanded"
		}
		logrus.Warn(detail)

		assessment.Violations = append(assessment.Violations, detail)
	}

	c.recordUpgradeEvent(assessment)
	return assessment, nil
}

// checkPVCMinSize determines whether a PVC was created from a
// VirtualMachineImage and, if so, whether its requested storage size is
// below that image's minimal disk size.
// It returns:
//   - (false, nil, nil) if PVC has no source image, or its size already
//     meets the minimum
//   - (true, minSize, nil) if PVC is undersized, along with the required
//     minimum size
//   - (false, nil, err) if the source image or its minimal disk size could
//     not be determined
func (c *Checker) checkPVCMinSize(pvc *corev1.PersistentVolumeClaim) (bool, *resource.Quantity, error) {
	image, err := util.GetPVCSourceImage(pvc, c.imageCache)
	if err != nil {
		return false, nil, err
	}
	if image == nil {
		return false, nil, nil
	}

	minSize, err := util.GetImageDiskSizeQuantity(image)
	if err != nil {
		return false, nil, err
	}

	if pvc.Spec.Resources.Requests.Storage().Cmp(*minSize) >= 0 {
		return false, nil, nil
	}

	return true, minSize, nil
}

func (c *Checker) recordUpgradeEvent(assessment *Assessment) {
	if c.upgradeName == "" {
		return
	}

	upgrade, err := c.upgradeCache.Get(util.HarvesterSystemNamespaceName, c.upgradeName)
	if err != nil {
		logrus.Warnf("failed to record image volume size check event: %v", err)
		return
	}

	eventType := corev1.EventTypeNormal
	message := fmt.Sprintf("Scanned %d volume(s), found %d smaller than their source image's virtual size.",
		assessment.Scanned, len(assessment.Violations))

	if len(assessment.Violations) > 0 {
		eventType = corev1.EventTypeWarning
		// Note, the event message has a length limitation, thus we can not
		// print the whole 'Violations' field here.
		message += " See upgrade-helper logs for details."
	}

	c.recorder.Event(upgrade, eventType, imageVolumeSizeCheckCompleted, message)
}
