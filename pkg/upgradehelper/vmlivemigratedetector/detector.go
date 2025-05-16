package vmlivemigratedetector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rancher/wrangler/v3/pkg/kv"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/retry"

	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"

	"github.com/harvester/harvester/pkg/util"
	"github.com/harvester/harvester/pkg/util/virtualmachineinstance"
)

type DetectorOptions struct {
	KubeConfigPath string
	KubeContext    string
	Shutdown       bool
	RestoreVM      bool
	NodeName       string
}

type VMLiveMigrateDetector struct {
	kubeConfig  string
	kubeContext string

	nodeName  string
	shutdown  bool
	restoreVM bool

	virtClient kubecli.KubevirtClient
}

func NewVMLiveMigrateDetector(options DetectorOptions) *VMLiveMigrateDetector {
	return &VMLiveMigrateDetector{
		kubeConfig:  options.KubeConfigPath,
		kubeContext: options.KubeContext,
		nodeName:    options.NodeName,
		shutdown:    options.Shutdown,
		restoreVM:   options.RestoreVM,
	}
}

func (d *VMLiveMigrateDetector) Init() (err error) {
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

	return
}

func (d *VMLiveMigrateDetector) Run(ctx context.Context) error {
	if d.nodeName == "" {
		return fmt.Errorf("please specify a node name")
	}

	// Get all VMs running on the specified node, except for the upgrade-related ones
	nodeReq, err := labels.NewRequirement("kubevirt.io/nodeName", selection.Equals, []string{d.nodeName})
	if err != nil {
		return err
	}
	notUpgradeReq, err := labels.NewRequirement("harvesterhci.io/upgrade", selection.DoesNotExist, nil)
	if err != nil {
		return err
	}
	selector := labels.NewSelector().Add(*nodeReq).Add(*notUpgradeReq)
	listOptions := metav1.ListOptions{
		LabelSelector: selector.String(),
	}
	vmiList, err := d.virtClient.VirtualMachineInstance("").List(ctx, listOptions)
	if err != nil {
		return err
	}
	vmis := make([]*kubevirtv1.VirtualMachineInstance, 0, len(vmiList.Items))
	for i := range vmiList.Items {
		vmis = append(vmis, &vmiList.Items[i])
	}

	// Get all nodes
	nodeList, err := d.virtClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	nodes := make([]*v1.Node, 0, nodeList.Size())
	for i := range nodeList.Items {
		nodes = append(nodes, &nodeList.Items[i])
	}

	nonLiveMigratableVMNames, err := virtualmachineinstance.GetAllNonLiveMigratableVMINames(vmis, nodes)
	if err != nil {
		return err
	}

	logrus.Infof("Non-migratable VM(s): %v", nonLiveMigratableVMNames)

	if d.shutdown {
		for _, namespacedName := range nonLiveMigratableVMNames {
			namespace, name := kv.RSplit(namespacedName, "/")
			if err := d.virtClient.VirtualMachine(namespace).Stop(ctx, name, &kubevirtv1.StopOptions{}); err != nil {
				return err
			}

			// if restoreVM is enabled, we need to add a label to the VM
			// to indicate that it should be restored after the upgrade
			if d.restoreVM {
				logrus.Infof("Adding restoreVM label to VM %s", namespacedName)

				payload, _ := json.Marshal([]util.PatchStringValue{
					{
						Op:    "add",
						Path:  "/metadata/labels/" + strings.ReplaceAll(util.LabelRestoreVMAfterUpgrade, "/", "~1"), // JSON Patch format requires '/' to be replaced with '~1'
						Value: d.nodeName,
					},
				})

				patchErr := retry.OnError(
					retry.DefaultBackoff,
					func(err error) bool { return err != nil && !apierrors.IsNotFound(err) },
					func() error {
						_, err := d.virtClient.VirtualMachine(namespace).Patch(ctx, name, types.JSONPatchType, payload, metav1.PatchOptions{})
						return err
					},
				)
				if patchErr != nil {
					logrus.Errorf("failed to patch VM %s with restoreVM label after retries: %v", namespacedName, patchErr)
					return patchErr
				}
			}

			logrus.Infof("vm %s was administratively stopped", namespacedName)
		}
	}

	return nil
}
