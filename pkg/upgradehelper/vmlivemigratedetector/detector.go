package vmlivemigratedetector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rancher/wrangler/v3/pkg/condition"
	"github.com/rancher/wrangler/v3/pkg/kv"
	"github.com/rancher/wrangler/v3/pkg/name"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
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
	harvNodeController "github.com/harvester/harvester/pkg/controller/master/node"
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
	RestoreVM      bool
	Upgrade        string
}

type VMLiveMigrateDetector struct {
	kubeConfig  string
	kubeContext string

	nodeName string
	shutdown bool

	restoreVM   bool
	upgradeName string

	virtClient   kubecli.KubevirtClient
	factory      *ctlharvester.Factory
	upgradeCache ctlharvesterv1.UpgradeCache
	recorder     record.EventRecorder
}

func NewVMLiveMigrateDetector(options DetectorOptions) *VMLiveMigrateDetector {
	return &VMLiveMigrateDetector{
		kubeConfig:  options.KubeConfigPath,
		kubeContext: options.KubeContext,
		nodeName:    options.NodeName,
		shutdown:    options.Shutdown,
		restoreVM:   options.RestoreVM,
		upgradeName: options.Upgrade,
	}
}

func (d *VMLiveMigrateDetector) Init() (err error) {
	if d.nodeName == "" {
		logrus.Fatal("please specify a node name")
	}
	if d.restoreVM && d.upgradeName == "" {
		logrus.Fatal("--upgrade {upgrade_name} must be specified when --restore-vm is used")
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

	factory, err := ctlharvester.NewFactoryFromConfig(restConfig)
	if err != nil {
		logrus.Fatalf("cannot obtain harvester factory: %v", err)
	}
	d.factory = factory
	d.upgradeCache = d.factory.Harvesterhci().V1beta1().Upgrade().Cache()

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

	if err := d.factory.Sync(ctx); err != nil {
		return fmt.Errorf("failed to sync factory: %w", err)
	}

	nodes, err := d.getAllNodes(ctx)
	if err != nil {
		return err
	}

	listOptions, err := d.getVMIListOptions(nodes)
	if err != nil {
		return err
	}

	vmis, err := d.getVMIs(ctx, listOptions)
	if err != nil {
		return err
	}

	nonLiveMigratableVMNames, err := getNonLiveMigratableVMNames(vmis, nodes)
	if err != nil {
		return err
	}

	logrus.Infof("Non-migratable VM(s): %v", nonLiveMigratableVMNames)

	if d.restoreVM {
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

func (d *VMLiveMigrateDetector) getAllNodes(ctx context.Context) ([]*v1.Node, error) {
	// filter out witness nodes as they are not available for vm migration
	nodeReq, err := labels.NewRequirement(harvNodeController.HarvesterWitnessNodeLabelKey, selection.DoesNotExist, nil)
	if err != nil {
		return nil, err
	}
	nodeList, err := d.virtClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: nodeReq.String()})
	if err != nil {
		return nil, err
	}
	nodes := make([]*v1.Node, 0, nodeList.Size())
	for i := range nodeList.Items {
		nodes = append(nodes, &nodeList.Items[i])
	}
	return nodes, nil
}

func (d *VMLiveMigrateDetector) getVMIs(ctx context.Context, opts metav1.ListOptions) ([]*kubevirtv1.VirtualMachineInstance, error) {
	vmiList, err := d.virtClient.VirtualMachineInstance("").List(ctx, opts)
	if err != nil {
		return nil, err
	}
	vmis := make([]*kubevirtv1.VirtualMachineInstance, 0, len(vmiList.Items))
	for i := range vmiList.Items {
		vmis = append(vmis, &vmiList.Items[i])
	}
	return vmis, nil
}

func (d *VMLiveMigrateDetector) getVMIListOptions(nodes []*v1.Node) (metav1.ListOptions, error) {
	// All VMs are non-migratable if there is only one node
	if len(nodes) == 1 {
		return metav1.ListOptions{}, nil
	}
	nodeReq, err := labels.NewRequirement("kubevirt.io/nodeName", selection.Equals, []string{d.nodeName})
	if err != nil {
		return metav1.ListOptions{}, err
	}
	notUpgradeReq, err := labels.NewRequirement("harvesterhci.io/upgrade", selection.DoesNotExist, nil)
	if err != nil {
		return metav1.ListOptions{}, err
	}
	return metav1.ListOptions{
		LabelSelector: labels.NewSelector().Add(*nodeReq).Add(*notUpgradeReq).String(),
	}, nil
}

func getNonLiveMigratableVMNames(vmis []*kubevirtv1.VirtualMachineInstance, nodes []*v1.Node) ([]string, error) {
	// All VMs are non-migratable if there is only one node
	if len(nodes) == 1 {
		names := make([]string, 0, len(vmis))
		for _, vmi := range vmis {
			names = append(names, vmi.Namespace+"/"+vmi.Name)
		}
		return names, nil
	}
	return virtualmachineinstance.GetAllNonLiveMigratableVMINames(vmis, nodes)
}

func getRestoreVMNames(vmis []*kubevirtv1.VirtualMachineInstance, nonMigratableVMNames []string) []string {
	restoreVMs := make([]string, 0, 0)

	// filter out paused VMs and upgrade repo VM from the non-migratable VMs
	excludeVMs := make(map[string]struct{})
	for _, vmi := range vmis {
		if vmPaused.IsTrue(vmi) || vmi.Labels["harvesterhci.io/upgrade"] != "" {
			namespacedName := fmt.Sprintf("%s/%s", vmi.Namespace, vmi.Name)
			excludeVMs[namespacedName] = struct{}{}
		}
	}
	for _, name := range nonMigratableVMNames {
		if _, exist := excludeVMs[name]; !exist {
			restoreVMs = append(restoreVMs, name)
		}
	}
	return restoreVMs
}
func (d *VMLiveMigrateDetector) getConfigMapName() string {
	return name.SafeConcatName(d.upgradeName, util.RestoreVMConfigMap)
}

func (d *VMLiveMigrateDetector) createOrUpdateConfigMap(ctx context.Context, restoreVMNames []string) error {
	vmNames := strings.Join(restoreVMNames, ",")
	name := d.getConfigMapName()
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
					newConfigMap := &v1.ConfigMap{
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
						fmt.Sprintf("ConfigMap %s/%s created", util.HarvesterSystemNamespaceName, d.getConfigMapName()))
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
