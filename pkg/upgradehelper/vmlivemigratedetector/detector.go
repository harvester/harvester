package vmlivemigratedetector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rancher/wrangler/v3/pkg/condition"
	"github.com/rancher/wrangler/v3/pkg/kv"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/retry"

	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"

	harvNodeController "github.com/harvester/harvester/pkg/controller/master/node"
	harvclient "github.com/harvester/harvester/pkg/generated/clientset/versioned"
	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
)

const (
	vmPaused condition.Cond = "Paused"
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

	nodeName  string
	shutdown  bool
	restoreVM bool
	upgrade   string

	virtClient kubecli.KubevirtClient
	harvClient harvclient.Interface
}

type PatchStringValue struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

func NewVMLiveMigrateDetector(options DetectorOptions) *VMLiveMigrateDetector {
	return &VMLiveMigrateDetector{
		kubeConfig:  options.KubeConfigPath,
		kubeContext: options.KubeContext,
		nodeName:    options.NodeName,
		shutdown:    options.Shutdown,
		restoreVM:   options.RestoreVM,
		upgrade:     options.Upgrade,
	}
}

func (d *VMLiveMigrateDetector) Init() (err error) {
	if d.restoreVM && d.upgrade == "" {
		logrus.Fatal("restoreVM is true while upgrade is not set")
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
		logrus.Fatalf("cannot obtain KubeVirt client: %v\n", err)
	}

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		logrus.Fatalf("cannot obtain rest config: %v\n", err)
	}

	d.harvClient, err = harvclient.NewForConfig(restConfig)
	if err != nil {
		logrus.Fatalf("cannot obtain harvester client: %v\n", err)
	}

	return
}

func (d *VMLiveMigrateDetector) Run(ctx context.Context) error {
	if d.nodeName == "" {
		return fmt.Errorf("please specify a node name")
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
		nodeUID := getNodeUID(d.nodeName, nodes)
		if nodeUID == "" {
			return fmt.Errorf("cannot find node %s UID", d.nodeName)
		}
		getRestoreVMNames := getRestoreVMNames(vmis, nonLiveMigratableVMNames)
		logrus.Infof("Patch upgrade CR with restoreVM annotation: %v", getRestoreVMNames)
		if err := d.patchUpgradeCR(ctx, getRestoreVMNames, nodeUID); err != nil {
			return err
		}
	}

	if d.shutdown {
		for _, namespacedName := range nonLiveMigratableVMNames {
			namespace, name := kv.RSplit(namespacedName, "/")
			if err := d.virtClient.VirtualMachine(namespace).Stop(ctx, name, &kubevirtv1.StopOptions{}); err != nil {
				return err
			}
			logrus.Infof("vm %s was administratively stopped", namespacedName)
		}
	}

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

func (d *VMLiveMigrateDetector) patchUpgradeCR(ctx context.Context, restoreVMNames []string, nodeUID string) error {
	annoKey := util.AnnotationRestoreVMPrefix + nodeUID
	annoValue := strings.Join(restoreVMNames, ",")
	payload, err := json.Marshal([]PatchStringValue{
		{
			Op: "add",
			// JSON Patch format requires '/' to be replaced with '~1'
			Path:  "/metadata/annotations/" + strings.ReplaceAll(annoKey, "/", "~1"),
			Value: annoValue,
		},
	})
	if err != nil {
		return err
	}

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		upgrade, err := d.harvClient.HarvesterhciV1beta1().Upgrades(util.HarvesterSystemNamespaceName).
			Get(ctx, d.upgrade, metav1.GetOptions{})
		if err != nil {
			return err
		}

		if value, ok := upgrade.Annotations[annoKey]; ok && value == annoValue {
			// already up-to-date
			return nil
		}

		_, err = d.harvClient.HarvesterhciV1beta1().Upgrades(util.HarvesterSystemNamespaceName).
			Patch(ctx, d.upgrade, types.JSONPatchType, payload, metav1.PatchOptions{})
		return err
	})
	return err
}

func getNodeUID(nodeName string, nodes []*v1.Node) string {
	for _, node := range nodes {
		if node.Name == nodeName {
			return string(node.UID)
		}
	}
	return ""
}
