package vmlivemigratedetector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rancher/wrangler/v3/pkg/condition"
	ctlcore "github.com/rancher/wrangler/v3/pkg/generated/controllers/core"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/kv"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"

	harvv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	ctlharvester "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
)

const (
	vmPaused condition.Cond = "Paused"

	RestoreVMConfigMapFailed  = "RestoreVMConfigMapFailed"
	RestoreVMConfigMapCreated = "RestoreVMConfigMapCreated"
	VMShutdownFailed          = "VMShutdownFailed"
	VMShutdownCompleted       = "VMShutdownCompleted"
)

type DetectorOptions struct {
	KubeConfigPath string
	KubeContext    string
	Shutdown       bool
	NodeName       string
	Upgrade        string
}

type VMLiveMigrateDetector struct {
	kubeConfig  string
	kubeContext string

	nodeName    string
	shutdown    bool
	upgradeName string

	virtClient   kubecli.KubevirtClient
	harvFactory  *ctlharvester.Factory
	upgradeCache ctlharvesterv1.UpgradeCache
	coreFactory  *ctlcore.Factory
	nodeCache    ctlcorev1.NodeCache
	recorder     record.EventRecorder
}

func NewVMLiveMigrateDetector(options DetectorOptions) *VMLiveMigrateDetector {
	return &VMLiveMigrateDetector{
		kubeConfig:  options.KubeConfigPath,
		kubeContext: options.KubeContext,
		nodeName:    options.NodeName,
		shutdown:    options.Shutdown,
		upgradeName: options.Upgrade,
	}
}

func (d *VMLiveMigrateDetector) Init() (err error) {
	if d.nodeName == "" {
		logrus.Fatal("please specify a node name")
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{
			ExplicitPath: d.kubeConfig,
		},
		&clientcmd.ConfigOverrides{
			ClusterInfo:    clientcmdapi.Cluster{},
			CurrentContext: d.kubeContext,
		},
	)

	d.virtClient, err = kubecli.GetKubevirtClientFromClientConfig(clientConfig)
	if err != nil {
		logrus.Fatalf("cannot obtain KubeVirt client: %v", err)
	}

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		logrus.Fatalf("cannot obtain rest config: %v", err)
	}

	harvFactory, err := ctlharvester.NewFactoryFromConfig(restConfig)
	if err != nil {
		logrus.Fatalf("cannot obtain harvester factory: %v", err)
	}
	d.harvFactory = harvFactory
	d.upgradeCache = harvFactory.Harvesterhci().V1beta1().Upgrade().Cache()

	coreFactory, err := ctlcore.NewFactoryFromConfig(restConfig)
	if err != nil {
		logrus.Fatalf("cannot obtain harvester factory: %v", err)
	}
	d.coreFactory = coreFactory
	d.nodeCache = coreFactory.Core().V1().Node().Cache()

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: d.virtClient.CoreV1().Events(util.HarvesterSystemNamespaceName)})
	d.recorder = broadcaster.NewRecorder(
		scheme.Scheme,
		corev1.EventSource{Component: "vm-live-migrate-detector", Host: d.nodeName},
	)

	return
}

func (d *VMLiveMigrateDetector) Run(ctx context.Context) error {
	defer func() {
		// wait for events to be flushed
		time.Sleep(10 * time.Second)
	}()

	if err := d.harvFactory.Sync(ctx); err != nil {
		return fmt.Errorf("failed to sync havester factory: %w", err)
	}
	if err := d.coreFactory.Sync(ctx); err != nil {
		return fmt.Errorf("failed to sync havester factory: %w", err)
	}

	vmis, err := d.getVMIs(ctx)
	if err != nil {
		return err
	}

	nodes, err := d.nodeCache.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	nonLiveMigratableVMNames, err := virtualmachineinstance.GetAllNonLiveMigratableVMINames(vmis, nodes)
	if err != nil {
		return err
	}

	logrus.Infof("Non-migratable VM(s): %v", nonLiveMigratableVMNames)

	if d.upgradeName != "" {
		vmNames := getRestoreVMNames(vmis, nonLiveMigratableVMNames)
		logrus.Infof("Store vm info to configmap: %v", vmNames)
		err = d.createOrUpdateConfigMap(ctx, vmNames)
		if err != nil {
			d.recordUpgradeEvent(corev1.EventTypeWarning, RestoreVMConfigMapFailed, err.Error())
			return err
		}
	}

	vmSuccessCnt := 0
	vmFailedCnt := 0
	if d.shutdown {
		for _, namespacedName := range nonLiveMigratableVMNames {
			namespace, name := kv.RSplit(namespacedName, "/")
			if err := d.virtClient.VirtualMachine(namespace).Stop(ctx, name, &kubevirtv1.StopOptions{}); err != nil {
				d.recordUpgradeEvent(corev1.EventTypeNormal, VMShutdownFailed,
					fmt.Sprintf("Shutdown failed for VM %s on node %s, error: %v", namespacedName, d.nodeName, err))
				logrus.Errorf("failed to stop VM %s: %v", namespacedName, err)
				vmFailedCnt++
			} else {
				vmSuccessCnt++
			}
			logrus.Infof("vm %s was administratively stopped", namespacedName)
		}
	}

	d.recordUpgradeEvent(corev1.EventTypeNormal, VMShutdownCompleted,
		fmt.Sprintf("Shutdown completed for %d VM(s) on node %s, success: %d, failed: %d ", len(nonLiveMigratableVMNames), d.nodeName, vmSuccessCnt, vmFailedCnt))

	return nil
}

func (d *VMLiveMigrateDetector) getVMIs(ctx context.Context) ([]*kubevirtv1.VirtualMachineInstance, error) {
	// filter out VMs that are not on the current node
	nodeReq, err := labels.NewRequirement("kubevirt.io/nodeName", selection.Equals, []string{d.nodeName})
	if err != nil {
		return nil, fmt.Errorf("failed to create node label requirement: %w", err)
	}
	options := metav1.ListOptions{LabelSelector: labels.NewSelector().Add(*nodeReq).String()}

	vmiList, err := d.virtClient.VirtualMachineInstance("").List(ctx, options)
	if err != nil {
		return nil, err
	}
	vmis := make([]*kubevirtv1.VirtualMachineInstance, 0, len(vmiList.Items))
	for i := range vmiList.Items {
		vmis = append(vmis, &vmiList.Items[i])
	}
	return vmis, nil
}

func getRestoreVMNames(vmis []*kubevirtv1.VirtualMachineInstance, candidateVMNames []string) []string {
	restoreVMs := make([]string, 0)

	// exclude paused VMs and upgrade repo VM from the candidate VMs
	// as they are not supposed to be restored
	excludeVMs := make(map[string]struct{})
	for _, vmi := range vmis {
		if vmPaused.IsTrue(vmi) || vmi.Labels["harvesterhci.io/upgrade"] != "" {
			namespacedName := fmt.Sprintf("%s/%s", vmi.Namespace, vmi.Name)
			excludeVMs[namespacedName] = struct{}{}
		}
	}
	for _, name := range candidateVMNames {
		if _, exist := excludeVMs[name]; !exist {
			restoreVMs = append(restoreVMs, name)
		}
	}
	return restoreVMs
}

func (d *VMLiveMigrateDetector) createOrUpdateConfigMap(ctx context.Context, restoreVMNames []string) error {
	vmNames := strings.Join(restoreVMNames, ",")
	name := util.GetRestoreVMConfigMapName(d.upgradeName)
	namespace := util.HarvesterSystemNamespaceName
	upgrade, err := d.upgradeCache.Get(util.HarvesterSystemNamespaceName, d.upgradeName)
	if err != nil {
		return fmt.Errorf("failed to get Upgrade CR %s: %w", d.upgradeName, err)
	}

	return retry.OnError(
		retry.DefaultBackoff,
		func(err error) bool {
			return errors.IsConflict(err) || errors.IsServerTimeout(err)
		},
		func() error {
			configMap, err := d.virtClient.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					// create a new ConfigMap if it does not exist
					newConfigMap := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									Name:       upgrade.Name,
									Kind:       "Upgrade",
									UID:        upgrade.UID,
									APIVersion: harvv1.SchemeGroupVersion.String(),
								},
							},
						},
						Data: map[string]string{
							d.nodeName: vmNames,
						},
					}
					_, createErr := d.virtClient.CoreV1().ConfigMaps(namespace).Create(ctx, newConfigMap, metav1.CreateOptions{})
					if createErr != nil && !errors.IsAlreadyExists(createErr) {
						return fmt.Errorf("failed to create ConfigMap: %w", createErr)
					}
					d.recordUpgradeEvent(corev1.EventTypeNormal, RestoreVMConfigMapCreated,
						fmt.Sprintf("ConfigMap %s/%s created", util.HarvesterSystemNamespaceName, util.GetRestoreVMConfigMapName(d.upgradeName)))
					return nil
				}
				return fmt.Errorf("failed to get ConfigMap: %w", err)
			}

			// update the existing ConfigMap
			if configMap.Data == nil {
				configMap.Data = map[string]string{}
			}
			configMap.Data[d.nodeName] = vmNames

			_, updateErr := d.virtClient.CoreV1().ConfigMaps(namespace).Update(ctx, configMap, metav1.UpdateOptions{})
			if updateErr != nil {
				return fmt.Errorf("failed to update ConfigMap: %w", updateErr)
			}
			return nil
		})
}

func (d *VMLiveMigrateDetector) recordUpgradeEvent(eventType, reason, message string) {
	if d.upgradeName == "" {
		return
	}
	upgrade, err := d.upgradeCache.Get(util.HarvesterSystemNamespaceName, d.upgradeName)
	if err != nil {
		logrus.Warnf("record event failed to get Upgrade CR %s: %v", d.upgradeName, err)
		return
	}
	logrus.Info("Recording event for upgrade", d.upgradeName, ":", eventType, reason, message)
	d.recorder.Event(upgrade, eventType, reason, message)
}
